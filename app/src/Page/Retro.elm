module Page.Retro
    exposing
        ( empty
        , mount
        , socketUpdate
        , update
        , view
        )

import Bulma
import Data.Column as Column
import Data.Retro as Retro
import Dict
import DragAndDrop
import EveryDict
import Html exposing (Html)
import Html.Attributes as Attr
import Page.RetroModel exposing (..)
import Page.RetroMsg exposing (Msg(..))
import Port
import Route
import Sock
import Views.Footer
import Views.Retro.Discussing
import Views.Retro.Header
import Views.Retro.Presenting
import Views.Retro.Thinking
import Views.Retro.Voting


empty : Model
empty =
    { retro = Retro.empty
    , inputs = EveryDict.empty
    , dnd = DragAndDrop.empty
    , lastRevealed = Nothing
    }


mount : String -> Sock.Sender Msg -> Cmd Msg
mount retroId sender =
    Sock.joinRetro sender retroId


update : Sock.Sender Msg -> Msg -> Model -> ( Model, Cmd Msg )
update sender msg model =
    case msg of
        Vote columnId cardId ->
            model ! [ Sock.vote sender columnId cardId ]

        Unvote columnId cardId ->
            model ! [ Sock.unvote sender columnId cardId ]

        SetStage stage ->
            { model | retro = Retro.setStage stage model.retro, lastRevealed = Nothing }
                ! [ Sock.stage sender (toString stage) ]

        Reveal columnId cardId ->
            { model | lastRevealed = Just cardId } ! [ Sock.reveal sender columnId cardId ]

        DnD subMsg ->
            case DragAndDrop.isDrop subMsg model.dnd of
                Just ( ( columnFrom, cardFrom ), ( columnTo, maybeCardTo ) ) ->
                    case model.retro.stage of
                        Retro.Thinking ->
                            if columnFrom /= columnTo then
                                { model | dnd = DragAndDrop.empty } ! [ Sock.move sender columnFrom columnTo cardFrom ]
                            else
                                model ! []

                        Retro.Voting ->
                            case maybeCardTo of
                                Just cardTo ->
                                    if cardFrom /= cardTo then
                                        { model | dnd = DragAndDrop.empty } ! [ Sock.group sender columnFrom cardFrom columnTo cardTo ]
                                    else
                                        model ! []

                                Nothing ->
                                    model ! []

                        _ ->
                            model ! []

                Nothing ->
                    { model | dnd = DragAndDrop.update subMsg model.dnd } ! []

        ChangeInput columnId input ->
            { model | inputs = EveryDict.insert columnId (String.trim input) model.inputs } ! []

        CreateCard columnId ->
            { model | inputs = EveryDict.remove columnId model.inputs }
                ! [ EveryDict.get columnId model.inputs
                        |> Maybe.map (Sock.add sender columnId)
                        |> Maybe.withDefault Cmd.none
                  ]

        DeleteCard columnId cardId ->
            model ! [ Sock.delete sender columnId cardId ]

        EditCard columnId cardId ->
            { model | retro = Retro.editingCard columnId cardId True model.retro } ! []

        UpdateCard columnId cardId contentId ->
            { model
                | inputs = EveryDict.remove columnId model.inputs
                , retro = Retro.editingCard columnId cardId False model.retro
            }
                ! [ Sock.edit sender contentId columnId cardId (EveryDict.get columnId model.inputs |> Maybe.withDefault "") ]

        DiscardEditCard columnId cardId ->
            { model | inputs = EveryDict.remove columnId model.inputs, retro = Retro.editingCard columnId cardId False model.retro } ! []

        Navigate route ->
            model ! [ Route.navigate route ]

        SignOut ->
            model ! [ Port.signOut () ]


parseStage : String -> Maybe Retro.Stage
parseStage s =
    case s of
        "Thinking" ->
            Just Retro.Thinking

        "Presenting" ->
            Just Retro.Presenting

        "Voting" ->
            Just Retro.Voting

        "Discussing" ->
            Just Retro.Discussing

        _ ->
            Nothing


socketUpdate : Maybe String -> Sock.Msg -> Model -> ( Model, Cmd Msg )
socketUpdate username msg model =
    case msg of
        Sock.Stage { stage } ->
            case parseStage stage of
                Just s ->
                    { model | retro = Retro.setStage s model.retro, lastRevealed = Nothing } ! []

                Nothing ->
                    model ! []

        Sock.Card { columnId, cardId, revealed, votes, totalVotes } ->
            let
                card =
                    { id = cardId
                    , votes = votes
                    , totalVotes = totalVotes
                    , revealed = revealed
                    , contents = []
                    , editing = False
                    }
            in
            { model | retro = Retro.addCard columnId card model.retro } ! []

        Sock.Content { contentId, columnId, cardId, author, cardText } ->
            let
                content =
                    { id = contentId
                    , text = cardText
                    , author = author
                    }
            in
            { model | retro = Retro.addContent columnId cardId content model.retro } ! []

        Sock.Column { columnId, columnName, columnOrder } ->
            let
                column =
                    { id = columnId, name = columnName, order = columnOrder, cards = EveryDict.empty }
            in
            { model | retro = Retro.addColumn column model.retro } ! []

        Sock.Move { columnFrom, columnTo, cardId } ->
            { model | retro = Retro.moveCard columnFrom columnTo cardId model.retro } ! []

        Sock.Reveal { columnId, cardId } ->
            { model | retro = Retro.revealCard columnId cardId model.retro, lastRevealed = Just cardId } ! []

        Sock.Group { columnFrom, cardFrom, columnTo, cardTo } ->
            { model | retro = Retro.groupCards ( columnFrom, cardFrom ) ( columnTo, cardTo ) model.retro } ! []

        Sock.Vote { userId, columnId, cardId } ->
            if Just userId == username then
                { model | retro = Retro.voteCard 1 columnId cardId model.retro } ! []
            else
                { model | retro = Retro.totalVoteCard 1 columnId cardId model.retro } ! []

        Sock.Unvote { userId, columnId, cardId } ->
            if Just userId == username then
                { model | retro = Retro.voteCard -1 columnId cardId model.retro } ! []
            else
                { model | retro = Retro.totalVoteCard -1 columnId cardId model.retro } ! []

        Sock.Delete { columnId, cardId } ->
            { model | retro = Retro.removeCard columnId cardId model.retro } ! []

        Sock.Error err ->
            Debug.log ("Sock.Error: " ++ toString err) model ! []

        _ ->
            model ! []


view : String -> Model -> Html Msg
view username model =
    Html.div [ Attr.class "site-content" ]
        [ Views.Retro.Header.view username model.retro.stage
        , Bulma.section [ Attr.class "fill-height x-auto-scroll" ]
            [ Html.div [ Attr.class "container is-fluid" ]
                [ case model.retro.stage of
                    Retro.Discussing ->
                        Views.Retro.Discussing.view model

                    Retro.Thinking ->
                        Views.Retro.Thinking.view username model

                    Retro.Presenting ->
                        Views.Retro.Presenting.view username model

                    Retro.Voting ->
                        Views.Retro.Voting.view model
                ]
            ]
        , Views.Footer.view
        ]
